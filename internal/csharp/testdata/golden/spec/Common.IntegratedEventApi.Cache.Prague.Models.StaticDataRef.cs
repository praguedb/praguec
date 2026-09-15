using MessagePack;

namespace Common.IntegratedEventApi.Cache.Prague.Models;

[MessagePackObject]
public class StaticDataRef
{
    [Key(0)]
    public long Id { get; set; }

    [Key(1)]
    public string Name { get; set; }

    [Key(2)]
    public string? ProviderSourceKey { get; set; }

    [Key(3)]
    public long ProviderKey { get; set; }
}
