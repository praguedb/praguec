using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(Prague.Models.BPTicketTypeCache))]
[DataCacheTopic("Cache.[v:brandId].TicketType")]
public sealed partial class TicketType
{
    [Key(0)]
    [DataCacheKey]
    public (long EventId, long TicketTypeId) Key => (EventId, TicketTypeId);

    [Key(1)]
    public long EventId { get; set; }

    [Key(2)]
    public long Id { get; set; }

    [IgnoreMember]
    public long TicketTypeId
    {
        get => Id;
        set => Id = value;
    }

    [Key(3)]
    public string Name { get; set; }

    [Key(6)]
    public TicketTypeStatus Status { get; set; }

    [Key(7)]
    public TicketTypeStatus? StatusManual { get; set; }

    [Key(8)]
    public TicketTypeStatus? StatusWatchDog { get; set; }

    [Key(21)]
    public decimal? LimitManualSingle { get; set; }

    [Key(24)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    [DataCacheFromTimestamp]
    public long Timestamp { get; set; }

    [Key(25)]
    public List<Quote> Quotes { get; set; }
}
