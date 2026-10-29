using Common.EventApi.Cache.Prague.Models;
using MessagePack;

namespace BrandReader.Services.Cache;

[MessagePackObject]
public sealed partial class SlimTicketType
{
    [Key(0)]
    [DataCacheKey]
    public (long EventId, long TicketTypeId) Id => (EventId, TicketTypeId);

    [Key(1)]
    public long EventId { get; set; }

    [Key(2)]
    public long TicketTypeId { get; set; }

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
    public long Timestamp { get; set; }

    [Key(25)]
    public List<SlimQuote> Quotes { get; set; }
}
