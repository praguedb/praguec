using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(Prague.Models.BPShowCache))]
[DataCacheTopic("Cache.[v:brandId].Show")]
public sealed partial class Show
{
    [Key(0)]
    [DataCacheKey]
    public long EventId { get; set; }

    [Key(1)]
    public long GenreId { get; set; }

    [Key(2)]
    public long CountryId { get; set; }

    [Key(3)]
    public long TourId { get; set; }

    [Key(4)]
    public long? HeadlinerId { get; set; }

    [Key(5)]
    public long? OpenerId { get; set; }

    [Key(6)]
    public Venue? Venue { get; set; }

    [Key(14)]
    public int? ListingId { get; set; }

    [Key(23)]
    public EventStatus EventStatus { get; set; }

    [Key(27)]
    public long EventDate { get; set; }

    [Key(34)]
    public long ProviderTimestamp { get; set; }

    [Key(35)]
    public decimal? LiveLimitManualSingle { get; set; }

    [Key(46)]
    public List<StaticDataRef> Performers { get; set; }

    [Key(54)]
    public StaticDataRef Genre { get; set; }

    [Key(55)]
    public StaticDataRef Country { get; set; }

    [Key(56)]
    public StaticDataRef Tour { get; set; }

    [Key(57)]
    public StaticDataRef? Headliner { get; set; }

    [Key(58)]
    public StaticDataRef? Opener { get; set; }
}
